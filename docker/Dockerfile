FROM alpine:3.20

RUN apk add --no-cache ca-certificates git

COPY knowing /usr/local/bin/knowing

ENTRYPOINT ["knowing"]
