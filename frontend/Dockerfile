# Build stage
FROM node:20-alpine AS builder

WORKDIR /app

# VITE_PHASE is baked into the JS bundle at build time by Vite.
# Pass it via docker-compose build args (see docker-compose.yml args section).
ARG VITE_PHASE=99
ENV VITE_PHASE=${VITE_PHASE}

COPY package.json ./
RUN npm install --legacy-peer-deps

COPY . .

RUN npm run build

# Runtime stage — nginx
FROM nginx:alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
