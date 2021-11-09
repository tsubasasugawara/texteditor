FROM golang:alpine

ENV ROOT=/go/src/app
WORKDIR ${ROOT}

ADD . .
RUN go mod init
