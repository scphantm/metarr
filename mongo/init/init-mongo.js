// Runs once, automatically, the first time the mongodb container starts
// with an empty data volume (see docker-entrypoint-initdb.d in the
// official mongo image docs). Provisions the "metarr" database with two
// application users:
//
//   - app       readWrite  -> user profiles + saved queries
//   - readonly  read       -> everything the Search screen touches
//
// The readonly user is the actual enforcement mechanism behind "no
// updates/deletes from the search screen": it physically cannot write,
// regardless of what the application sends it.

const dbName = process.env.MONGO_DATABASE || "metarr";
const appUser = process.env.MONGO_APP_USERNAME || "app";
const appPassword = process.env.MONGO_APP_PASSWORD || "apppassword";
const readonlyUser = process.env.MONGO_READONLY_USERNAME || "readonly";
const readonlyPassword = process.env.MONGO_READONLY_PASSWORD || "readonlypassword";

const targetDb = db.getSiblingDB(dbName);

targetDb.createUser({
  user: appUser,
  pwd: appPassword,
  roles: [{ role: "readWrite", db: dbName }],
});

targetDb.createUser({
  user: readonlyUser,
  pwd: readonlyPassword,
  roles: [{ role: "read", db: dbName }],
});

// Seed an empty media collection so the readonly user has something to
// query against out of the box.
targetDb.createCollection("media");
