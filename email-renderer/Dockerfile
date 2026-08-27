FROM mcr.microsoft.com/playwright:v1.62.0-noble

WORKDIR /app

COPY package.json package-lock.json ./
RUN PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm ci --omit=dev --no-audit --no-fund

# Keep every local ES module alongside the entry point. This prevents a
# successful Node syntax test on the host from becoming a missing-module crash
# only after the production image starts.
COPY *.mjs ./

USER pwuser

EXPOSE 8080

CMD ["node", "server.mjs"]
