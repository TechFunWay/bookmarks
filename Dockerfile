FROM scratch

WORKDIR /app

ARG TARGETARCH
ARG VERSION
COPY release/${VERSION}/bookmarks-${VERSION}-linux-${TARGETARCH}/bookmarks /app/bookmarks
COPY release/${VERSION}/bookmarks-${VERSION}-linux-${TARGETARCH}/reset-password /app/reset-password

EXPOSE 8901

VOLUME /app/data

CMD ["/app/bookmarks", "-deviceType=docker"]
