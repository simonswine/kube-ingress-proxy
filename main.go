package main

import (
	log "github.com/Sirupsen/logrus"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/util/intstr"
)

const appName = "kube-inress-proxy"
const appVersion = "0.0.1"

func main() {

	log.Infof("Starting of %s %s", appName, appVersion)

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
