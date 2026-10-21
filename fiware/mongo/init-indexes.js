// Entity indexes recommended by Orion's performance-tuning guide (doc/manuals/admin/perf_tuning.md).
// Orion creates no indexes on the entities collection itself (only 2dsphere/TTL when needed).
// With -multiservice, entities of fiware-service <S> live in database "orion-<S>".
const service = process.env.FIWARE_SERVICE || "openiot";
const dbNames = ["orion", "orion-" + service];

for (const name of dbNames) {
  const ents = db.getSiblingDB(name).entities;
  ents.createIndex({ "_id.servicePath": 1, "_id.id": 1, "_id.type": 1 });
  ents.createIndex({ creDate: 1 });
  ents.createIndex({ "_id.id": 1 });
  ents.createIndex({ "_id.type": 1 });
  ents.createIndex({ "_id.servicePath": 1 });
  ents.createIndex({ attrNames: 1 });
  print(`indexes ensured on ${name}.entities`);
}
