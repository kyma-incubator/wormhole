FROM scratch

ADD wormhole-dispatcher /wormhole-dispatcher
ADD dispatcher.pem /dispatcher.pem
ADD dispatcher-key.pem /dispatcher-key.pem

ENTRYPOINT ["/wormhole-dispatcher"]
