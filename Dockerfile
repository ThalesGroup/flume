FROM golang:1.11.1-alpine

# ENV http_proxy=$http_proxy
# ENV http_proxy=http://$HTTP_PROXY
RUN apk --no-cache add make git curl bash fish

# build tools
COPY ./Makefile /gosrc/
WORKDIR /gosrc
RUN make tools

COPY ./ /gosrc

CMD make all

# unset proxy vars, just used in build
# ENV http_proxy=
