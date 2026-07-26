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

## Future Roadmap

These are features I want to put into the system.

### Versioned save

When documents are saved they can be saved with cache time to live or permanent versioned typed

If the cached_ttl field is set it just sits there.  A scheduled job scans the documents and deletes
any documents from the database that exceed the ttl.

Versioned save is different.
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
I want to have a read/write agent that can be run on remote machines.  This agent can connect to the server via the Redis caches

But the idea is that reading drives over the network is I/O expensive.  If I had a small as possible agent that could be run
on the physical nas machine, it can do all the file read/write operations locally and simply send the results over the event system
instead of reading the file system over the network, like NFS or whatever.  

In theory, this should give a big boost in speed to read/write operations.

The agent can be used for download operations as well, basically the equivalent of running wget from your machine over 
a network drive or locally on that server.  I'm thinking this for operations like image downloads.  That agent will 
be written in Go or Rust because we want them tiny and as fast as possible.  These languages are binary languages and are far faster than interpreted
languages like python, Java, C#, etc.

https://towardsdev.com/file-writing-speed-battle-node-python-go-rust-php-and-c-8a1c35ad870e
