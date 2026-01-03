FROM golang:1.25-alpine

ENV AIR_VERSION=v1.63.0 \
    GOLANGCI_LINT_VERSION=v2.7.2 \
    MIGRATE_VERSION=v4.19.1 \
    MOCKERY_VERSION=v3.6.1

RUN apk add --no-cache \
    git \
    make \
    curl \
    postgresql-client \
    tzdata \
    libc6-compat \
    netcat-openbsd

    
RUN go install github.com/air-verse/air@${AIR_VERSION}

RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@${MIGRATE_VERSION}

RUN go install github.com/vektra/mockery/v3@${MOCKERY_VERSION}

RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
    sh -s -- -b /go/bin v${GOLANGCI_LINT_VERSION}

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY ./docker/init.sh /usr/local/bin/init.sh
RUN chmod +x /usr/local/bin/init.sh

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]