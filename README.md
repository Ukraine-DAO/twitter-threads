# Twitter thread archiver/renderer

This piece of code is designed to run periodically, fetch new tweets from
configured threads and update generated Markdown files for each threads.

## How to use

All threads are listed in [`config.yml`](config.yml) under `root` key:

```yaml
root:
    my_thread: 123456789123456789
```

Key under `root` will be the name of the page (`my_thread.md`) and the value is
the ID of a tweet that belongs to the desired thread.

You can also specify a title for a thread:

```yaml
root:
    my_thread:
        thread_id: 123456789123456789
        title: My fancy thread
```

To group threads into a tree structure you can use `subdirs`:

```yaml
root:
    my_thread: 123456789123456789
    subdirs:
        important:
            my_important_thread: 987654321987654321
```

Keys on the first level under `subdirs` will become directories, and under each
of them you can specify threads just like under `root` (in fact, `root` is
actually defined as a subdir, so it behaves identically).

Definitive information on all knobs available is in
[`common/config.go`](common/config.go).

### Which tweet ID to use?

If you put the ID of the first tweet in a thread, collector will try to fetch
the full tree of replies by the thread author and pick the longest chain out of
it.

If you put ID of a tweet further down the thread, collector will walk up from it
to the start of the thread *ignoring any branches*, and attempt to fetch the
full tree below the specified tweet.

Due to limitations of Twitter API, for older threads putting ID of the first
tweet might not work. But putting the ID of the last tweet should always work.

### Running locally

To run the generator locally you'll need:

1. Working Go compiler
2. Any valid Twitter bearer token (obtainable by
registering and creating an app on Twitter developer portal), stored in
`.secrets/twitter_bearer_token` file.
3. GNU `make`

First, update the generated files to their current state:

```sh
git submodule update --init --remote
```

Then run the collector and renderer:

```sh
make collect render
```

If it doesn't spit out anything looking like an error, you should get the output
in `generated/` directory.

## Technical details

For our purposes a thread is defined as the longest chain of replies by the same
user that starts from a tweet by that user.

To track that, collector stores the whole tree of replies by the thread author
in `state` branch.

Additionally, if the specified tweet ID is not the first in the thread, branches
that start off from tweets before it are ignored, i.e., we fetch only a single
chain of tweets there by following `replied_to` relationship.

### Limitations

Without Academic Research access tweets search endpoint only returns tweets from
the last 7 days, which isn't very useful.

So instead of that, we're fetching tweets specified in the config by ID (which
works regardless of tweet age), and then fetch the timeline of the tweet author.
Timeline returns up to 3200 last tweets, which usually covers a lot more than
7 days.

Reconstructing threads from user's timeline is not particularly hard given that
`conversation_id` is the same for all tweets in a thread and `replied_to` points
to the previous tweet.

To avoid fetching all 3200 tweets every time, we store the point up to which we
fetched the timeline the last time in `state` branch, so the next time we can
fetch only new tweets after it. Whenever a new thread by the same user is added
that value gets reset in order to have a chance to fetch tweets belonging to the
newly added thread.

