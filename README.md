## github-artifact-fetcher

Github Artifact Fetcher is a web app written in Go reacting to Github Actions Webhooks and fetching artifacts produced by the received Action run.

This project supports [distributhor/workflow-webhook@v2](https://github.com/marketplace/actions/workflow-webhook-action) action.
Take a look at example configuration for your webhook step

```yaml
- name: fetcher webhook
  uses: distributhor/workflow-webhook@v2
  env:
    webhook_url: http://example.com/receiver
    webhook_secret: My$up3Rp@s$w0rD
    webhook_type: json
    data: '{"run_id": ${{ github.run_id }}}'
```

Make sure that `run_id` is defined like in the example above as it's crucial for the fetcher.

#### How does it work?

Fetcher starts a web server on a corresponding host:port (see Configuration section) and reacts to exactly one endpoint which is also configurable (default: `/receive`).

When your action reaches the Webhook step it sends a payload to fetcher, e.g.

```json
{
  "event":"push",
  "repository":"vatsimnerd/simwatch",
  "commit":"93cc5f9ed2844237c6c169a3ca3761a726a4b930",
  "ref":"refs/heads/gh-actions",
  "head":"",
  "workflow":"Simwatch",
  "data":{
    "run_id":2431090188
  },
  "requestID":"72efa85b-811d-4c05-8101-2c715bcd43a8"
}
```

Fetcher checks if the `repository` and `workflow` fields match fetcher artifacts configuration and if it does, makes a request to Github API to find URLs for the artifacts of the current `run_id`, downloads zip archive and unzips it to the destination folder configured for this artifact. That's it.

There are lifecycle callbacks for each artifact downloaded, `before` and `after` which may be configured for each artifact independently. Some useful variables are available for these callbacks:

```
GITHUB_ARTIFACT_ID
GITHUB_ARTIFACT_NAME
GITHUB_ARTIFACT_SIZE
GITHUB_ARTIFACT_URL
GITHUB_ARTIFACT_DOWNLOAD_URL
```

#### Configuration

Configuration is quite self-explanatory

```yaml
addr: localhost:5000
endpoint: /receive
artifacts:
  - name: simwatch-server
    repo: vatsimnerd/simwatch
    path: "."
    github_token: xxxxSECRETxxxx
    filter:
      event: push
      workflow: simwatch
    after:
      - echo Name is $GITHUB_ARTIFACT_NAME
```
