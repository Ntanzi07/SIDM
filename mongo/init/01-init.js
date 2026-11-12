// Executado pelo Mongo apenas na PRIMEIRA vez que o volume é criado.
db = db.getSiblingDB("sidm");

db.createCollection("images");      // TODO: adicionar validador $jsonSchema
db.createCollection("image_history");
db.createCollection("codes");

db.images.createIndex({ equipment: 1, captured_at: -1 });
db.images.createIndex({ camera: 1, captured_at: -1 });
db.images.createIndex({ day_time: 1, captured_at: -1 });
db.images.createIndex({ weather: 1, captured_at: -1 });

db.image_history.createIndex({ image_id: 1, changed_at: -1 });
