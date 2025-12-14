# This is a renovate-friendly source of Docker images.
FROM python:3.13.5-slim-bullseye@sha256:5b9fc0d8ef79cfb5f300e61cb516e0c668067bbf77646762c38c94107e230dbc AS python
FROM otel/weaver:v0.20.0@sha256:fa4f1c6954ecea78ab1a4e865bd6f5b4aaba80c1896f9f4a11e2c361d04e197e AS weaver
FROM avtodev/markdown-lint:v1@sha256:6aeedc2f49138ce7a1cd0adffc1b1c0321b841dc2102408967d9301c031949ee AS markdown
