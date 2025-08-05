FROM node:22-alpine

WORKDIR /app

# Build-time environment variables
ARG VITE_API_URL
ARG VITE_ALLOWED_REDIRECT_DOMAINS
ARG VITE_ALLOWED_HOSTS
ARG VITE_DEFAULT_DOMAINS

# Set them as environment variables for build
ENV VITE_API_URL=$VITE_API_URL
ENV VITE_ALLOWED_REDIRECT_DOMAINS=$VITE_ALLOWED_REDIRECT_DOMAINS
ENV VITE_ALLOWED_HOSTS=$VITE_ALLOWED_HOSTS
ENV VITE_DEFAULT_DOMAINS=$VITE_DEFAULT_DOMAINS

# Copy and install dependencies
COPY package*.json ./
RUN npm install

# Kaynak kodu kopyala
COPY . .

# Build the application
RUN npm run build

# Port
EXPOSE 80

# Set API_URL with environment variable
ENV API_URL=http://api:3000/api/v1

# Serve static files with Vite - remove all host restrictions
CMD ["npm", "run", "preview", "--", "--host", "0.0.0.0", "--port", "80", "--strictPort", "false"] 