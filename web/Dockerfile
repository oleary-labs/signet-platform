FROM node:20-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci

FROM node:20-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# NEXT_PUBLIC_* values are inlined at build time, so they are build args rather
# than runtime environment. Every one the app reads needs a line here — one
# that is only set at runtime reaches the browser as undefined, which fails as
# a feature that silently does nothing rather than as an error.
ARG NEXT_PUBLIC_API_URL=http://localhost:8088
ARG NEXT_PUBLIC_SITE_URL=http://localhost:3000
ARG NEXT_PUBLIC_GOOGLE_CLIENT_ID=
ARG NEXT_PUBLIC_BUNDLER_URL=
ARG NEXT_PUBLIC_USE_PAYMASTER=false
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_SITE_URL=$NEXT_PUBLIC_SITE_URL
ENV NEXT_PUBLIC_GOOGLE_CLIENT_ID=$NEXT_PUBLIC_GOOGLE_CLIENT_ID
ENV NEXT_PUBLIC_BUNDLER_URL=$NEXT_PUBLIC_BUNDLER_URL
ENV NEXT_PUBLIC_USE_PAYMASTER=$NEXT_PUBLIC_USE_PAYMASTER
RUN npm run build

FROM node:20-alpine
WORKDIR /app
ENV NODE_ENV=production
RUN addgroup -g 10001 signet && adduser -D -u 10001 -G signet signet
# .next, not .next-build: `npm run build` writes to the canonical directory and
# `npm start` reads from it. Setting NEXT_DIST_DIR in either would break this
# copy, and the failure is a build-time "no source files" with no hint why.
COPY --from=build /app/.next ./.next
COPY --from=build /app/public ./public
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/package.json ./package.json
USER signet
EXPOSE 3000
CMD ["npm", "run", "start"]
