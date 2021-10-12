FROM golang:1.16.7
LABEL PROJECT="kconfig-deployer"

COPY src /go/src/kconfig-deployer/
WORKDIR /go/src/kconfig-deployer/

RUN go get -d

RUN go install

RUN mkdir -p bin
RUN go build -o bin/

ENTRYPOINT ["/go/src/konfig-deployer/bin/kconfig-deployer"]