# syntax=docker/dockerfile:1
FROM golang:1.16.4-stretch
RUN go get -u github.com/beego/bee
ENV GO111MODULE=on
ENV GOFLAGS=-mod=vendor
WORKDIR app
EXPOSE 8010
CMD ["bee", "run"]