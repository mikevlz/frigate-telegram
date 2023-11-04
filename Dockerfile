FROM golang:1.21.3-alpine3.18 AS builder

WORKDIR $GOPATH/frigate-telegram
COPY . $GOPATH/frigate-telegram

RUN apk --no-cache add binutils
RUN go build .
RUN strip frigate-telegram

FROM alpine:3.18

RUN apk --no-cache add tzdata

COPY --from=builder /go/frigate-telegram/frigate-telegram /frigate-telegram

USER 1000

ENTRYPOINT ["/frigate-telegram"]
