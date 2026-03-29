FROM busybox:1.37
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
RUN mkdir -p /workspace /tmp && chown 1000:1000 /workspace /tmp
USER 1000:1000
ENTRYPOINT ["/bin/sh", "/entrypoint.sh"]
