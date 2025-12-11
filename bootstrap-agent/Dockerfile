FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /src/bootstrap-agent

FROM docker:26-cli

WORKDIR /opt/citizen

COPY --from=build /src/bootstrap-agent /usr/local/bin/bootstrap-agent

RUN apk add --no-cache git

ENV BOOTSTRAP_BIND="0.0.0.0:8085" \
    BOOTSTRAP_DATA_DIR="/opt/citizen/data"

EXPOSE 8085

ENTRYPOINT ["/usr/local/bin/bootstrap-agent"]
