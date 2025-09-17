# Build and serve the React application
FROM node:18-alpine

# Set working directory
WORKDIR /app

# Copy package.json and package-lock.json
COPY package*.json ./
COPY vite.config.js ./
COPY tailwind.config.js ./
COPY postcss.config.js ./

# Install dependencies
RUN npm install

# Copy the rest of the application code
COPY . .

# Accept build-time arguments
ARG VITE_API_BASE
ENV VITE_API_BASE=$VITE_API_BASE

# Build the application
RUN npm run build

# Expose port 80
EXPOSE 80

# Start the server
CMD ["npm", "start"]