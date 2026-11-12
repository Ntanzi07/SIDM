// Executado pelo Mongo apenas na PRIMEIRA vez que o volume é criado.
db = db.getSiblingDB("sidm");

db.createCollection("images");     // { _id: sha1, package, status: pending|verified, ... }
db.createCollection("image_history");

// Pacotes: cada imagem pertence a UM pacote (pasta dataset/<nome>/).
db.createCollection("packages");    // { name, created_by, created_at }
db.packages.createIndex({ name: 1 }, { unique: true });

db.createCollection("cameras");     // { name: "hikvision-233", active: true }  (name já sanitizado)
db.createCollection("equipments");  // { name: "8003-MF4739", active: true }    (name já sanitizado)

db.cameras.createIndex({ name: 1 }, { unique: true });
db.equipments.createIndex({ name: 1 }, { unique: true });

db.images.createIndex({ package: 1, status: 1, queued_at: 1 });  // fila de trabalho e progresso de cada pacote
db.images.createIndex({ equipment: 1, captured_at: -1 });
db.images.createIndex({ camera: 1, captured_at: -1 });
db.images.createIndex({ day_time: 1, captured_at: -1 });
db.images.createIndex({ weather: 1, captured_at: -1 });

db.image_history.createIndex({ image_id: 1, changed_at: -1 });
