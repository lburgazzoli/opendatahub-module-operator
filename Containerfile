# Build all real module operator binaries and package them into one image.
FROM registry.access.redhat.com/ubi10/go-toolset:latest AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=0.0.0-dev
ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
ARG GIT_REPO=unknown
ARG EXCLUDED_MODULES="opendatahub-mymodule-operator"

USER 0
WORKDIR /workspace

# Copy the full repo because the root image discovers modules dynamically.
COPY . .

RUN bash hack/scripts/build-all-modules.sh && chmod 0755 hack/scripts/run-module.sh

# Use UBI 10 minimal so the launcher can stay a small POSIX shell script.
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest
WORKDIR /
COPY --from=builder /out/bin/ /opt/odh-modules/bin/
COPY --from=builder /out/manifests/ /opt/odh-modules/manifests/
COPY --from=builder /out/modules.txt /opt/odh-modules/modules.txt
COPY --from=builder /workspace/hack/scripts/run-module.sh /usr/local/bin/run-module
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/run-module"]
