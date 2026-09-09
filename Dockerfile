FROM    golang:1.26-alpine AS builder
RUN     apk add --no-cache make git
WORKDIR /app
COPY    . ./
RUN     make build

FROM        alpine:3.22 AS production
ENV         USER_UID="1000" \
            USER_GID="1000" \
            USER_NAME="mittwald-container-action"

RUN         apk add --no-cache ca-certificates && \
            addgroup -g ${USER_GID} ${USER_NAME} && \
            adduser -D -u ${USER_UID} -G "${USER_NAME}" "${USER_NAME}"

COPY        --from=builder /app/mittwald-container-action /mittwald-container-action

USER        ${USER_NAME}

ENTRYPOINT  ["/mittwald-container-action"]
