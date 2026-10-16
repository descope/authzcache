# Load-test image only. Built from a locally cross-compiled binary (see build.sh) so it needs
# no GitHub token. Alpine rather than scratch so `docker exec` works for RSS inspection.
FROM alpine:3

COPY app /app

EXPOSE 8189 6060
ENTRYPOINT ["/app"]
