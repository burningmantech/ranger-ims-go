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

## TestUsers

When working locally, you might want to configure a simple set of IMS users,
so you don't need a full Clubhouse DB. To do this, copy `conf/testusers.example.go`
to `conf/testusers.go` (that'll make it actually load as the server starts).
In that file, you'll configure the dummy users you want for your Directory.

After that, you just need to toggle on the TestUsers feature in your `.env`:

```shell
IMS_DIRECTORY="TestUsers"
```
