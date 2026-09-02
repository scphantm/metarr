---
name: implement-design
description: Implements a design from the Matt Pocock workflow
disable-model-invocation: true
---

# Setup

Make sure the session is connected to a PR. If there isn't a currently assigned PR, stop and ask the user to create one.

All output from this will be tuned for token efficiency.

## Steps

The issue being passed will be the design issue. Assign that issue to the current PR so its closed when the PR is
closed.

The issue will have sub issues under it. loop thru each of those sub issues one at a time running the `/implement` skill
in a new clear session.

When they are completed, assign the issue to the PR for auto close and move on to the next issue until complete.
