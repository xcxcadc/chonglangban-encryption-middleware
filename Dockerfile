FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/chonglangban-middleware .

FROM alpine:3.20
RUN addgroup -S middleware && adduser -S middleware -G middleware
WORKDIR /app
COPY --from=build /out/chonglangban-middleware /app/chonglangban-middleware
COPY .env.example /app/.env.example
USER middleware
EXPOSE 3000
ENTRYPOINT ["/app/chonglangban-middleware"]
