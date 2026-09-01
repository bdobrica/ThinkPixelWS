# Service container image

The root `Dockerfile` builds the `thinkpixelws` service as a statically linked,
trimmed Go binary and copies only that binary into a distroless runtime. Both
the official Go builder and distroless runtime are pinned by multi-platform OCI
digest. The runtime has no shell or package manager and runs as UID/GID 65532.

Build it with:

```sh
make image
```

The safe process defaults bind to loopback. A container deployment must opt in
to externally reachable listeners, for example:

```sh
docker run --rm --read-only --cap-drop=ALL \
  -e THINKPIXELWS_HTTP_LISTEN_ADDRESS=0.0.0.0:8080 \
  -e THINKPIXELWS_METRICS_LISTEN_ADDRESS=0.0.0.0:9090 \
  -p 127.0.0.1:8080:8080 -p 127.0.0.1:9090:9090 \
  thinkpixelws:dev
```

The service writes logs to standard output and needs no writable filesystem for
the current baseline. Production orchestration should keep the root filesystem
read-only, drop all Linux capabilities, prevent privilege escalation, and place
the metrics listener behind deployment network policy. Secrets remain external
references and must not be copied into the image or supplied as build arguments.

`make check-service-image` enforces digest pins, static compilation, the numeric
non-root user, and the exec-form entrypoint. Image vulnerability scanning, SBOM,
signature, and provenance remain release-packaging gates tracked later in the
release plan.
