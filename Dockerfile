FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cineroom ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/cineroom /app/cineroom
COPY web /app/web
ENV ADDR=:8080 DATA_DIR=/data APP_ORIGIN=http://localhost:8080 MAX_UPLOAD_MB=1024
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/app/cineroom"]
