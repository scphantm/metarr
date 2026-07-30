# Code Quality
* All code should have descriptive variable names and follow standard accepted clean code practices.

# API Changes
On any changes made to the api, reflect those same changes on the cli application as well

# Changes to the configuration file structure

Any time changes are made to the configuration file structure, also execute these commands

* update the config api to include crud methods for the new configuration settings in the configuration router
* update the database initialization in `/cmd/api/main.go` to initialize the setting.