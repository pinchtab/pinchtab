# Custom fixtures image: nginx + baked-in test config.
#
# We bake the nginx config into the image instead of bind-mounting it,
# so the e2e suite is immune to "did docker actually pick up the new
# volume" failure modes when iterating locally or in CI.
#
# Static fixture *content* (HTML/JS/etc.) is still bind-mounted at
# /usr/share/nginx/html so it can be edited without rebuilding.
FROM nginx:alpine

# Replace the stock default site with our test-aware config that adds
# httpbin-compatible /redirect/N, /redirect-to, and /get endpoints.
COPY default.conf /etc/nginx/conf.d/default.conf

# Build-time guarantees:
# 1. config syntactically valid (catches typos)
# 2. our new endpoints are actually present (catches "wrong file copied")
# 3. no second default_server is hiding in the image elsewhere (catches
#    a stale image with leftover stock config taking precedence)
RUN nginx -t \
 && grep -q 'location = /redirect-to'          /etc/nginx/conf.d/default.conf \
 && grep -q 'location = /redirect-to-internal' /etc/nginx/conf.d/default.conf \
 && grep -q 'location = /redirect/1'           /etc/nginx/conf.d/default.conf \
 && grep -q 'location = /get'                  /etc/nginx/conf.d/default.conf
