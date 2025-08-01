# IMS Config

The IMSConfig struct (see imsconfig.go) holds all the configuration
for an IMS server. The settings are loaded into this struct through
the following path:

1. Start with the values returned by `conf.DefaultIMS()`
2. Override that with any values in `${RepoRoot}/.env`
3. Override that with environment variables

The `.env` part is optional, but we assume that local development
will primarily make use of a `.env` file. There's an example version
already in the repo, so just do the following, then tweak the resultant
file to meet your needs.

```shell
cd "${git rev-parse --show-toplevel}"
cp .env-example .env
```
