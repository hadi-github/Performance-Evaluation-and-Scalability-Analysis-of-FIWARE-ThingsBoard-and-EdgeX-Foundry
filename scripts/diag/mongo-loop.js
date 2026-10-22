// sidecar (mongo image, mongosh): opcounters every 5 s as JSON lines. With PROFILE_RATE set: once the update rate
// has been >= PROFILE_RATE for three samples, wait 30 s, profile all application DBs (level 2) for 60 s, then print
// the aggregation by (ns, op, planSummary) and every collection's indexes.
const rate = Number(process.env.PROFILE_RATE || 0);
const appDbs = () => db.adminCommand({ listDatabases: 1 }).databases.map((d) => d.name)
  .filter((n) => !['admin', 'local', 'config'].includes(n));
let prev = null, okRuns = 0, profiled = false;
while (true) {
  const op = db.serverStatus().opcounters;
  const t = Date.now();
  print(JSON.stringify({ t_ms: t, opcounters: op }));
  if (rate > 0 && !profiled && prev) {
    const up = (op.update - prev.op.update) / ((t - prev.t) / 1000);
    okRuns = up >= rate ? okRuns + 1 : 0;
    if (okRuns >= 3) {
      sleep(30000);
      const dbs = appDbs();
      for (const n of dbs) {
        const d = db.getSiblingDB(n);
        d.setProfilingLevel(0); d.system.profile.drop();
        d.createCollection('system.profile', { capped: true, size: 256 * 1024 * 1024 });
      }
      const on = Date.now();
      for (const n of dbs) db.getSiblingDB(n).setProfilingLevel(2);
      sleep(60000);
      for (const n of dbs) db.getSiblingDB(n).setProfilingLevel(0);
      const off = Date.now();
      const agg = [];
      for (const n of dbs) {
        db.getSiblingDB(n).system.profile.aggregate([
          { $group: { _id: { ns: '$ns', op: '$op', plan: '$planSummary' }, n: { $sum: 1 }, millis: { $sum: '$millis' },
                      docsExamined: { $sum: '$docsExamined' }, keysExamined: { $sum: '$keysExamined' },
                      nreturned: { $sum: '$nreturned' }, nModified: { $sum: '$nModified' } } }]).forEach((r) => agg.push(r));
      }
      const idx = {};
      for (const n of dbs) {
        const d = db.getSiblingDB(n);
        for (const c of d.getCollectionNames().filter((c) => !c.startsWith('system.'))) idx[n + '.' + c] = d[c].getIndexes();
      }
      print(JSON.stringify({ profile: { on_ms: on, off_ms: off, dbs: dbs, groups: agg, indexes: idx } }));
      profiled = true;
    }
  }
  prev = { op: op, t: t };
  sleep(5000);
}
