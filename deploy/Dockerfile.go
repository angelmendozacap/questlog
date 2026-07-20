# Builds any backend/cmd/* binary. CMD_PATH selects which one (public-api | admin-api | migrate).
FROM golang:1.26-alpine AS build
ARG CMD_PATH
WORKDIR /src
COPY go.work go.work
COPY backend/go.mod backend/go.sum backend/
COPY backend/tools/go.mod backend/tools/
RUN cd backend && go mod download
COPY backend/ backend/
RUN cd backend && CGO_ENABLED=0 go build -o /out/app ./${CMD_PATH}

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
