# 普段の開発に使うツールをまとめた Dev Container。ツールの版は CI (.github/workflows) と magefiles/magefile.go に合わせる。
# ゲームの Mod を組み立てるときだけ使うもの (.NET SDK・MinGW・Python など) は入れず、mage package:<game> がその都度コンテナで動かす。
FROM golangci/golangci-lint:v2.15.0 AS golangci
FROM koalaman/shellcheck:v0.11.0 AS shellcheck
FROM rhysd/actionlint:1.7.12 AS actionlint
FROM zricethezav/gitleaks:v8.30.1 AS gitleaks

FROM mcr.microsoft.com/devcontainers/go:2-1.27-trixie
COPY --from=golangci /usr/bin/golangci-lint /usr/local/bin/golangci-lint
COPY --from=shellcheck /bin/shellcheck /usr/local/bin/shellcheck
COPY --from=actionlint /usr/local/bin/actionlint /usr/local/bin/actionlint
COPY --from=gitleaks /usr/bin/gitleaks /usr/local/bin/gitleaks
RUN GOBIN=/usr/local/bin GOFLAGS=-mod=mod GOPATH=/tmp/gopath GOCACHE=/tmp/gocache \
      go install github.com/magefile/mage@v1.17.2 \
    && rm -rf /tmp/gopath /tmp/gocache
