# Minimal Go build for the expense-api stand-in. Single static binary,
# distroless runtime, in-memory state.
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/expense-api ./...

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/expense-api /usr/local/bin/expense-api
EXPOSE 8093
ENTRYPOINT ["/usr/local/bin/expense-api"]
