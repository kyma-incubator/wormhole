FROM alpine:latest

LABEL name=wormhole-connector

# set UID=1000, GID=1000 to avoid running with root
ARG UNAME=wc
ARG UID=1000
RUN adduser -g $UNAME -u $UID -D $UNAME
USER $UNAME

# wormhole-connector should have a writable work directory.
WORKDIR /tmp

# We need to have entrypoint.sh included in the image, to be able to
# have a correct local address being passed to wormhole-connector.
ADD aci/entrypoint.sh /entrypoint.sh
ADD wormhole-connector /wormhole-connector

# Expose necessary ports to make it accessible by external containers:
# 1111 for Serf, 1112 for Raft, and 8080 for HTTPS.
EXPOSE 1111 1112 8080

ENTRYPOINT /entrypoint.sh
