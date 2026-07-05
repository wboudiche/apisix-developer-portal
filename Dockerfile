# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /portal ./cmd/portal

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /portal /portal
EXPOSE 8080
ENTRYPOINT ["/portal"]
