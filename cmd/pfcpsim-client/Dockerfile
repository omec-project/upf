# Stage mock-smf-build: builds the mockSMF docker image
FROM golang AS mock-smf-build
WORKDIR /mock-smf

COPY go.mod ./go.mod
COPY go.sum ./go.sum

RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 go build -o /bin/mock-smf cmd/pfcpsim-client/main.go

# Stage mock-smf: runtime image of mockSMF
FROM alpine AS mock-smf
COPY --from=mock-smf-build /bin/mock-smf /bin
ENTRYPOINT [ "/bin/mock-smf", "-r", "127.0.0.1", "-v", "-i",  "eth0" ]