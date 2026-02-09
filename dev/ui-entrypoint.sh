#!/bin/sh
set -e

# Replace the default API URL with the configured one in all JS bundles
if [ -n "$API_BASE_URL" ] && [ "$API_BASE_URL" != "http://localhost:8150" ]; then
  echo "Replacing API base URL with: $API_BASE_URL"
  find /usr/share/nginx/html/assets -name "*.js" -type f -exec \
    sed -i "s|http://localhost:8150|${API_BASE_URL}|g" {} +
fi

# Execute the original entrypoint
exec /docker-entrypoint.sh "$@"
