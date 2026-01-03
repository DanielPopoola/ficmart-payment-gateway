FROM golang:1.25-alpine

ENV AIR_VERSION=v1.63.0 \
    GOLANGCI_LINT_VERSION=v2.7.2 \
    MIGRATE_VERSION=v4.19.1 \
    MOCKERY_VERSION=v3.6.1

RUN apk add --no-cache git postgresql-client tzdata curl libc6-compat

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

RUN go install github.com/air-verse/air@${AIR_VERSION}

RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin ${GOLANGCI_LINT_VERSION}


RUN curl -L https://github.com/golang-migrate/migrate/releases/download/${MIGRATE_VERSION}/migrate.linux-amd64.tar.gz | tar xvz && \
    mv migrate /usr/local/bin/migrate

RUN go install github.com/vektra/mockery/v3@${MOCKERY_VERSION}


COPY ./docker/init.sh /usr/local/bin/init.sh
RUN chmod +x /usr/local/bin/init.sh

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]