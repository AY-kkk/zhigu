# syntax=docker/dockerfile:1
FROM node:22 AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ARG VITE_INTEL_ENABLED=true
ENV VITE_INTEL_ENABLED=$VITE_INTEL_ENABLED
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.27-alpine
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
ARG VERSION=dev
LABEL org.opencontainers.image.revision=${VCS_REF} \
      org.opencontainers.image.source=zhigu \
      org.opencontainers.image.created=${BUILD_DATE} \
      org.opencontainers.image.version=${VERSION}
USER 101
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 8080 8443
