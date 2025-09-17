# Build and serve the React application
FROM node:18-alpine as builder

# Set working directory
WORKDIR /app

# Copy files
COPY . ./

# Install dependencies
RUN npm install

<<<<<<< HEAD
# Copy the rest of the application code
COPY . .

# Accept build-time arguments
ARG VITE_API_BASE
ENV VITE_API_BASE=$VITE_API_BASE

=======
>>>>>>> f1790372dcb9ce58a72e00068b8899692c57fd7c
# Build the application
RUN npm run build && ls -la

FROM nginx:1.29-alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY tools/nginx.template.conf /etc/nginx/nginx.template.conf

RUN chmod -R 777 /etc/nginx /var/cache/nginx /var/run
EXPOSE 27182

USER nginx

# Start the server
CMD /bin/sh -c "envsubst '\
    \$STATE_MANAGER_URL' \
    < /etc/nginx/nginx.template.conf \
    > /etc/nginx/conf.d/default.conf" \
    && nginx -g 'daemon off;'