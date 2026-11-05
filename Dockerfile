FROM    golang:1.26.10-alpine3.24@sha256:a2012b4a1d1f9f7e9f44e0d39b805a901e856b1dd80b4e6ee4338db547050308 AS builder
RUN     apk add --no-cache make git
WORKDIR /app
COPY    . ./
RUN     make build

FROM        alpine:3.22.7@sha256:2e0b81d60597f449baa56dc25d355367869d2dca4cd57e866f1e2b52af3eb55f AS production
ENV         USER_UID="1000" \
            USER_GID="1000" \
            USER_NAME="mittwald-container-action"

RUN         apk add --no-cache ca-certificates && \
            addgroup -g ${USER_GID} ${USER_NAME} && \
            adduser -D -u ${USER_UID} -G "${USER_NAME}" "${USER_NAME}"

COPY        --from=builder /app/mittwald-container-action /mittwald-container-action

USER        ${USER_NAME}

ENTRYPOINT  ["/mittwald-container-action"]
