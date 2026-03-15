FROM scratch

WORKDIR /app

ARG TARGETARCH
COPY release/bookmarks-v1.9.0-linux-${TARGETARCH}/bookmarks /app/bookmarks

EXPOSE 8901

VOLUME /app/data

CMD ["/app/bookmarks"]