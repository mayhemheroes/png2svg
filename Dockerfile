FROM golang:1.19.1-buster as builder

RUN DEBIAN_FRONTEND=noninteractive \
	apt-get update && apt-get install -y build-essential 

ADD . /png2svg
WORKDIR /png2svg
RUN go mod tidy
WORKDIR /png2svg/cmd/png2svg
RUN go build

FROM golang:1.19.1-buster
COPY --from=builder /png2svg/cmd/png2svg/png2svg /
COPY --from=builder /png2svg/testdata /testsuite/

ENTRYPOINT ["/png2svg", "@@"]
