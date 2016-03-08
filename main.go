package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/util/intstr"
)

type IngressProxy struct {
	Ingress   *extensions.Ingress
	HttpPort  int16
	HttpsPort int16
	backends  map[string]*httputil.ReverseProxy
}

func NewIngressProxy() *IngressProxy {
	i := &IngressProxy{
		HttpPort:  8080,
		HttpsPort: 8443,
	}
	i.backends = make(map[string]*httputil.ReverseProxy)
	return i
}

func (ip *IngressProxy) urlFromBackend(b *extensions.IngressBackend) *url.URL {
	return &url.URL{
		Host: fmt.Sprintf(
			"%s.svc.%s.cluster.local:%d",
			b.ServiceName,
			ip.Ingress.ObjectMeta.Namespace,
			b.ServicePort.IntVal,
		),
		Scheme: "http",
	}
}

func (ip *IngressProxy) routeRequest(r *http.Request) *httputil.ReverseProxy {

	backend := ip.routeRequestToBackend(r)
	backendKey := fmt.Sprintf("%s:%s", backend.ServiceName, backend.ServicePort)

	if backendProxy, ok := ip.backends[backendKey]; ok {
		return backendProxy
	}

	backendProxy := httputil.NewSingleHostReverseProxy(
		ip.urlFromBackend(backend),
	)
	ip.backends[backendKey] = backendProxy

	return backendProxy
}

func (ip *IngressProxy) routeRequestToBackend(r *http.Request) *extensions.IngressBackend {

	for _, rule := range ip.Ingress.Spec.Rules {
		if strings.ToLower(rule.Host) != strings.ToLower(r.Host) {
			//skip if hostname does not match
			continue
		}

		matchingLen := 0
		var matchingBackend *extensions.IngressBackend
		matchingBackend = nil
		for _, path := range rule.HTTP.Paths {
			if strings.HasPrefix(r.URL.Path, path.Path) && len(path.Path) > matchingLen {
				matchingLen = len(path.Path)
				matchingBackend = &path.Backend
			}
		}
		if matchingBackend != nil {
			return matchingBackend
		}

	}

	return ip.Ingress.Spec.Backend
}

func (ip *IngressProxy) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-KubeIngressProxy", "go alter!")
	log.Infof("host=%s path=%s method=%s", r.Host, r.URL.Path, r.Method)
	ip.routeRequest(r).ServeHTTP(w, r)
}

func (ip *IngressProxy) Start() {
	http.HandleFunc("/", ip.handle)
	log.Infof("Start listening for HTTP on port %d", ip.HttpPort)
	http.ListenAndServe(fmt.Sprintf(":%d", ip.HttpPort), nil)
}

func main() {
	config := &extensions.Ingress{
		ObjectMeta: api.ObjectMeta{
			Name:      "ingress1",
			Namespace: "default",
		},
		Spec: extensions.IngressSpec{
			Backend: &extensions.IngressBackend{
				ServiceName: "service1",
				ServicePort: intstr.FromInt(8080),
			},
			Rules: []extensions.IngressRule{
				extensions.IngressRule{
					Host: "www.test.de",
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								extensions.HTTPIngressPath{
									Path: "/",
									Backend: extensions.IngressBackend{
										ServiceName: "service2",
										ServicePort: intstr.FromInt(8080),
									},
								},
							},
						},
					},
				},
				extensions.IngressRule{
					Host: "www.test.co.uk",
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								extensions.HTTPIngressPath{
									Path: "/",
									Backend: extensions.IngressBackend{
										ServiceName: "service3",
										ServicePort: intstr.FromInt(8080),
									},
								},
								extensions.HTTPIngressPath{
									Path: "/backend",
									Backend: extensions.IngressBackend{
										ServiceName: "service4",
										ServicePort: intstr.FromInt(8080),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ip := NewIngressProxy()
	ip.Ingress = config

	ip.Start()
}
