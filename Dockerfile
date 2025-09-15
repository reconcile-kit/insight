# Build and serve the React application
FROM node:18-alpine as builder

# Set working directory
WORKDIR /app

# Copy files
COPY . ./

# Install dependencies
RUN npm install

# Build the application
RUN npm run build && ls -la && tree

FROM nginx:1.29-alpine

COPY --from=builder /app/build/bundle /usr/share/nginx/html
COPY tools/nginx.template.conf /etc/nginx/nginx.template.conf

RUN chmod -R 777 /etc/nginx /var/cache/nginx /var/run
EXPOSE 80

USER nginx

# Start the server
CMD /bin/sh -c "envsubst '\
    \$STATE_MANAGER_URL' \
    < /etc/nginx/nginx.template.conf \
    > /etc/nginx/conf.d/default.conf" \
    && nginx -g 'daemon off;'