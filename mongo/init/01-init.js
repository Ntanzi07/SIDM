// Executado pelo Mongo apenas na PRIMEIRA vez que o volume é criado.
db = db.getSiblingDB("sidm");

db.createCollection("images");
db.createCollection("image_history");

db.createCollection("cameras");     // { name: "hikvision-233", active: true }  (name já sanitizado)
db.createCollection("equipments");  // { name: "8003-MF4739", active: true }    (name já sanitizado)

db.cameras.createIndex({ name: 1 }, { unique: true });
db.equipments.createIndex({ name: 1 }, { unique: true });

db.images.createIndex({ equipment: 1, captured_at: -1 });
db.images.createIndex({ camera: 1, captured_at: -1 });
db.images.createIndex({ day_time: 1, captured_at: -1 });
db.images.createIndex({ weather: 1, captured_at: -1 });

db.image_history.createIndex({ image_id: 1, changed_at: -1 });
