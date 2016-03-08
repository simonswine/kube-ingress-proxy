package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"k8s.io/kubernetes/pkg/apis/extensions"
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
		matchingBackend := -1

		for pos, path := range rule.HTTP.Paths {
			fullPath := r.URL.Path
			prefixPath := path.Path
			if strings.HasPrefix(fullPath, prefixPath) && len(prefixPath) > matchingLen {
				matchingLen = len(path.Path)
				matchingBackend = pos
			}
		}

		if matchingBackend != -1 {
			return &rule.HTTP.Paths[matchingBackend].Backend
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
