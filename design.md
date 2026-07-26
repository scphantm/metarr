# Overview

create the scafolding of the application with these parameters

* The base language for this application will be typescript for the front end and python for the backend.
* build an initial UI using https://github.com/Sportarr/Sportarr as the starting point.
* The server side will be written in python with a rest api done in FastAPI at https://pypi.org/project/fastapi/.
* Build a docker file that packages the entire application into a single docker file using the latest alpine node image
* create a docker compose file that creates an instance of MongoDB
* put the connection information for the backend services to connect to the database in a configuration file at the root of the project

## Search
The backend is in MongoDB.  The search page will allow you to enter mongodb queries and search your media library.

* Build a system that allows you to save your queries to your user profile in the database
* The user interface will be split in half.  The top half will be a query entry box
* The bottom screen will display the results of the query.  
* Very strict controls will be in place to prevent updates and deletes from this screen.  This will be query only.

Build the scaffold for a web api with the following requirements
* The language is the latest version of GoLang
* The primary data store is MongoDB
* Redis is used for the pub/sub, caching, and event driven backend systems

The system architecture will have both a pub/sub queue using Redis as the message queue and
an event driven backend using Redis as the backend server.  

all messages that flow in the backend will have correlation id's

There will be a heartbeat api.  This heartbeat will register an event on the queue.  A listener on that event queue will reply with the current date time and the correlation id. 
This is the response sent to the web client.  This is a blocking call

The second api will trigger a long running job.  The api will have the following requirements
* post to `api/tasks/sonarr_cache_data` with the body `{"command": "run"}` 
* The service will fire the event `sonarr_cache_data` on the event bus in a non-blocking way.
* The api will return to the client once the event is fired.
* On the backend, the SonarrCacheData listener sees the event, fires, and logs to the log file "event fired"


on the backend, build out a system capable of running long running jobs.  This system will mantain a queue of jobs to be run.  Develop a 
class structure in python to make creating new jobs easier.  To create a job, you extend the class and fill in the code for the run_job method. 
A rest api jobs/queue will take a string parameter.  That string parameter will map to a dictionary in the service, that will use the key passed 
to queue the correct job in the long running system.

Migrate the health check to run via the event queue.  the job queue convert to a redis pub sub queue to handle long running jobs


Add an application config system with the following requirements.
* The app config structure's is exampled in app_config.local.yaml.  build a model for this file so it can be stored in the mongo database.
* The model and rest of the code should be built with the assumption that there will only ever be one copy of this config document in the database
* Add an `api/config` get to the route that will retrieve the app config from mongo and return it to the web client.
* a put to `api/config` should update the configuration
* when the put comes in.  The service will fire an event `system_config_update` with the updated document as the payload and that will be picked up by a listener.  
    That listener will update the record in the mongo database.  Then it will update the global singleton configuration value to the new settings.