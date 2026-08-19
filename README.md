# Metarr

In our media collections, the media files themselves are obviously important.  But as your collection grows,
what becomes ever increasingly important is the maintenance of the metadata of your collection.  Its this metadata
that ensures things get named right in systems, play correctly, and sort and organize your collection.  

All the downloaders of the ServArr suite support writing metadata.  But that capability is limited by the number of 
sources they use and different formats.  Stack more systems on top of your media library like TV Tuners, image systems
and the like, and maintaining the metadata becomes nearly impossible.  TinyMediaManager does an ok, job, but it is very
slow and doesn't read tags from Sonarr or Radarr.  

Metarr is designed to maintain the metadata of your media collection so it can ultimatly be consumed
by Jellyfin.  (Other outputs possible).  

Unlike other *arr's, the database in this project is more for caching data from external systems than
metadata storage.  With Metarr, the final system of record for your metadata are the nfo files in the file system.  

## What this does not do

This does not look for missing media in your collection.  Other systems are designed to do that.  

## Local URI's

* [Swagger API](http://localhost:8080/swagger/index.html)
* [Mongo Express](http://10.0.0.22:6969/)
* [Redis Insights](http://localhost:5540/)

## Architecture

Metarr is two binaries.

**metarr-server** owns the API, MongoDB, and orchestration. It never touches the
media library — it has no filesystem access to it at all.

**metarr-agent** is a small static binary deployed next to the storage, on the
NAS itself where possible. It does every filesystem operation: walking
libraries, reading NFO files, inspecting artwork. It connects to Redis and
nothing else, holds no database credentials, and cannot open a database
connection — a test walks the build graph on every run to keep that true.

An agent is configured locally with two things: how to reach Redis, and its own
name. Everything else is published to it over Redis by the server. Start one and
it appears under **System > Agents** as connected but unconfigured; configuring
it means mapping the libraries the server knows about onto the paths that
machine knows them by. Records are always stored under the server's paths, so
the library reads the same however many agents produced it.

Agents run on Windows, Linux or macOS. Build one for a target with
`make dist`, which emits static binaries for each.

```
make run-server                                    # the API
make run-agent METARR_AGENT_SLUG=nas-01            # an agent alongside it
docker compose up                                  # both, plus Mongo and Redis
```

## Architectural features
Extensive use of caching.  I/O operations are the slowest operations in all of computer science.  And this application
does thousands of them.  To prevent you from rate limiting your API keys, and to cut back on compilation times, this system
caches virtually everything.  All the time to lives are configurable in the configuration system

Event driven backend.  Because I/O is so time expensive, Metarr takes advantage of an Event Driven Backend to spread jobs across
several cores in parallel.  This means many things happen at once, instead of
sequentially like other systems you may be used to.  Metarr follows a model called Eventually Consistent.  Meaning when you
hit save, the change you make may not be instantaneous.  Many things happen in parallel in the background and all have to finish
before the write actually happens.  Not when you click the save button.  Eventually Consistent means it will get there and update
just maybe not instantly

## UI

The UI will consist of 3 critical pieces.  
* Searches
* Workflows
* Automations

Searches will use [React Query Builder](https://react-querybuilder.js.org/docs/utils/import#custom-operators) to query the mongodb. 

From there, you will be able to build out workflows using [React Workflow](https://reactflow.dev/ui/components/animated-svg-edge) that are basically what i 
currently do with claude scripts

Automations are mapping search results, workflows, and some kind of trigger.  cron, event bus hook, webhook, etc and what agent to run on

Things like what i just wrote as a downloader will become workflows.  Thats going to be my first big task, taking my youtube downloader and making
it a workflow in metarr.

Workflows will be dry-run only to begin with.  There will be some kind of combination lock that puts the system into write mode that will
allow workflows being worked on to write.

metadata management itself will include a scraper system.  the one that grabs from all the different areas.

## List manager
Want a system that lets you compare my database with mdblists to group things, see what i already have, retag, request missing, etc.

Lists could be an input to workflows.  That way you can build workflows that process lists.

## Future Roadmap

These are features I want to put into the system.

### Versioned save

When documents are saved they can be saved with cache time to live or permanent versioned typed

If the cached_ttl field is set it just sits there.  A scheduled job scans the documents and deletes
any documents from the database that exceed the ttl.
ioned save is different.
* Save the initial document
* On update, it does a compare with the existing document and records the difference somehow.
* the get will have an overload, if you request a previous version, the service will use the history to reconstruct
    that old version of the file

### Safe rename
I want a system where you can run reports on different naming conventions to see what would change.  When you come up
with a standard pattern for naming your media, it should do the rename, AND update the correct darr to the new path
so things line up.  It should update jellyfin too if the api has the ability.

I want the rename system to have conditionals.  Like scripts.  Maybe Lua?  I don't know.  But the rename will not be 
a simple regex pattern, it will have conditionals.

### git style diff
show a diff between what metarr believes the it should change like git

### moment in time snapshot
I want to snapshot the metadata library.  That way i can compare what changed over time.

### Backup
I want to backup the metadata library.  all of it, everything except the video files themselves.

### Agents

Done — see Architecture above. The remaining work is to move download operations
(artwork fetching, and the wget-equivalent for external assets) onto the agent
as well, so those also happen next to the storage rather than across the network.

### Metadata proxy.
everything here is cached and versioned.  I want to build this so all other systems can point to it and
they get cached results and versioned so you can see how a document changed over a period of time.  That 
data will be used along side our data just like any other.

### Metadata update
This should have a way to update the other metadata registries if its possible thru their api.

### Poster manager
Think Kometa poster manager, you can build one or more poster and image profiles for a media item.  those poster bundles
will be stored somewhere and swapped in at will.  The different poster places will be huge for that.

# References
This system interfaces other systems.  These were the specifications that were used in building Metarr

## Directory Structures
* https://jellyfin.org/docs/general/server/media/shows/
* https://jellyfin.org/docs/general/server/media/movies

## Naming Conventions
* https://support.plex.tv/articles/naming-and-organizing-your-movie-media-files/
* https://support.plex.tv/articles/naming-and-organizing-your-tv-show-files/

## NFO Format
https://kodi.wiki/view/NFO_files/Templates