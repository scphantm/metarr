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
//
// It also seeds the singleton application config document (see
// internal/appconfig/model.go) with a freshly generated API key for each
// access-level category. Without this, there's no way to obtain the very
// first admin key needed to call PUT /api/config, since that route itself
// requires an admin key.

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

const appConfigCollection = targetDb.getCollection("app_config");

if (appConfigCollection.countDocuments({ _id: "app_config" }) === 0) {
  const adminKey = crypto.randomUUID();
  const userKey = crypto.randomUUID();
  const webhookKey = crypto.randomUUID();
  const readOnlyKey = crypto.randomUUID();

  appConfigCollection.insertOne({
    _id: "app_config",
    api_keys: {
      admin: [{ name: "Administrator Key", api_key: adminKey }],
      user: [{ name: "User Key", api_key: userKey }],
      webhook: [{ name: "Webhook Key", api_key: webhookKey }],
      read_only: [{ name: "Read Only Key", api_key: readOnlyKey }],
    },
    interfaces: { sonarr: [] },
  });

  print("");
  print("==================================================================");
  print("Metarr: generated default API keys (shown only once, save these):");
  print("  admin:     " + adminKey);
  print("  user:      " + userKey);
  print("  webhook:   " + webhookKey);
  print("  read_only: " + readOnlyKey);
  print("==================================================================");
  print("");
} else {
  print("Metarr: app_config document already exists, skipping API key seed.");
}
