# Build the React app, then serve the static bundle with nginx.
FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
# Template is rendered at startup via envsubst (API_UPSTREAM), then nginx serves
# the SPA and proxies /api to the apiserver.
COPY default.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
ENV API_UPSTREAM=http://wordpress-apiserver.wordpress-system.svc.cluster.local:80
EXPOSE 80
