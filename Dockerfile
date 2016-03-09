FROM golang:1.5

RUN go get github.com/tools/godep

WORKDIR /go/src/github.com/simonswine/kube-ingress-proxy/
ADD *.go ./
ADD Godeps ./Godeps/

RUN godep go test && godep go build

CMD ./kube-ingress-proxy
