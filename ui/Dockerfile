FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npx nx build admin-ui --configuration=production

FROM nginx:alpine3.21
RUN rm /etc/nginx/conf.d/default.conf && \
    chown -R nginx:nginx /var/cache/nginx /var/run /run
COPY --chown=nginx:nginx --from=build /app/dist/admin-ui/browser /usr/share/nginx/html
COPY --chown=nginx:nginx nginx.conf /etc/nginx/conf.d/default.conf
USER nginx
EXPOSE 80
