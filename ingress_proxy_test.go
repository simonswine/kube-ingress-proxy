package main

import (
	"testing"
	"net/http"
	"net/url"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/util/intstr"
)

func TestSampleConfigRouting(t *testing.T) {
	i := exampleIngress()

	r := http.Request{}
	r.Host = "www.test.de"
	r.URL = &url.URL{Path: "/any/page/asd"}
	b := i.routeRequestToBackend(&r)
	if b.ServiceName != "service2" {
		t.Errorf("request=%+v routed to wrong backend=%+v", r, b)
	}

	r = http.Request{}
	r.Host = "www.test.co.uk"
	r.URL = &url.URL{Path: "/any/page/asd"}
	b = i.routeRequestToBackend(&r)
	if b.ServiceName != "service3" {
		t.Errorf("request=%+v routed to wrong backend=%+v", r, b)
	}

	r = http.Request{}
	r.Host = "www.test.co.uk"
	r.URL = &url.URL{Path: "/backend/asd"}
	b = i.routeRequestToBackend(&r)
	if b.ServiceName != "service4" {
		t.Errorf("request=%+v routed to wrong backend=%+v", r, b)
	}

}

func exampleIngress() *IngressProxy {

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

	return ip
}
