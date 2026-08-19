# Planning

Claude is not permitted to edit any code under any circumstances without presenting a plan and having it approved first.  No exceptions.

# Code Quality
* All code should have descriptive variable names and follow standard accepted clean code practices.

# Changes to the configuration file structure

Any time changes are made to the configuration file structure, also execute these commands

* update the config api to include crud methods for the new configuration settings in the configuration router
* update the database initialization in `/cmd/api/main.go` to initialize the setting.

# Data update methods

When adding crud methods to the api for a block of data, instead of creating separate POST and PUT methods, 
create a single POST that upserts

# External API Calls

All http calls to external api's should be done with the cached http client.  This ensures that all calls are cached
and reusable based on the configuration settings.