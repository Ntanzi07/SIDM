# --- build ---
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /sidm ./cmd/server

# --- runtime ---
FROM alpine:3
COPY --from=build /sidm /sidm
EXPOSE 8080
ENTRYPOINT ["/sidm"]
