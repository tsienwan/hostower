# syntax=docker/dockerfile:1
# 多阶段构建：产物是一个静态二进制，放进没有 shell、没有包管理器的镜像里，非 root 运行。

FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
# 自我更新的来源和发布公钥：发布流程传进来；不传就是一个不会自我更新的构建。
ARG UPDATE_REPO="" UPDATE_KEY=""
# PREBUILT=1：直接用 dist/hostower_linux_<架构>（发布流程已经编译好、加了构建封签的那份），不在这里重新编译。
ARG PREBUILT=""
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
# 先编译增强采集程序（两种架构都要：Hostower 可能管着和自己架构不同的主机），它会被编译进 Hostower 里。
RUN mkdir -p /out/data && if [ "$PREBUILT" = 1 ]; then \
      install -m 755 "dist/hostower_linux_$TARGETARCH" /out/hostower; \
    else \
      go run ./tools/buildagent "$VERSION" && \
      CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
      go build -trimpath -ldflags "-s -w -X main.version=$VERSION -X main.updateRepo=$UPDATE_REPO -X main.updateKey=$UPDATE_KEY" -o /out/hostower ./cmd/hostower; \
    fi

# 宿主机版（docker build --target host）：容器部署也能开箱管理所在的这台机器。
# 它要以特权容器运行并进入宿主机的命名空间，等于宿主机上的 root；用法和取舍见 deploy/compose.host.yaml。
FROM alpine:3.22 AS host
# 自带一份静态的 docker 命令：宿主机上只有 Docker 服务、没有命令行时用它（宿主机自己有的优先）。
COPY --from=docker:29-cli /usr/local/bin/docker /opt/hostower/bin/docker
COPY --from=build /out/hostower /hostower
COPY --from=build /out/data /data
ENV HOSTOWER_DATA=/data \
    HOSTOWER_CONFIG=/config \
    HOSTOWER_LISTEN=0.0.0.0:18443 \
    HOSTOWER_LOCAL=host
EXPOSE 18443
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/hostower", "healthcheck"]
ENTRYPOINT ["/hostower"]
CMD ["serve"]

# 默认镜像（最后一个阶段）：不带 shell，非 root，管不到宿主机。
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hostower /hostower
COPY --from=build --chown=65532:65532 /out/data /data
ENV HOSTOWER_DATA=/data \
    HOSTOWER_CONFIG=/config \
    HOSTOWER_LISTEN=0.0.0.0:18443
USER 65532:65532
EXPOSE 18443
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/hostower", "healthcheck"]
ENTRYPOINT ["/hostower"]
CMD ["serve"]
