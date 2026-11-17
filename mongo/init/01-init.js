// Executado pelo Mongo apenas na PRIMEIRA vez que o volume é criado.
db = db.getSiblingDB("sidm");

db.createCollection("images");     // { _id: sha1, package, status: pending|verified, ... }
db.createCollection("image_history");

// Pacotes: cada imagem pertence a UM pacote (pasta dataset/<nome>/).
db.createCollection("packages");    // { name, created_by, created_at }
db.packages.createIndex({ name: 1 }, { unique: true });

// Perfis de import: o "modelo" de cada formato de CSV ({ name, spec: "<json>", ... }).
db.createCollection("import_profiles");
db.import_profiles.createIndex({ name: 1 }, { unique: true });

db.createCollection("cameras");     // { name: "hikvision-233", active: true }  (name já sanitizado)
db.createCollection("equipments");  // { name: "8003-MF4739", active: true }    (name já sanitizado)

db.cameras.createIndex({ name: 1 }, { unique: true });
db.equipments.createIndex({ name: 1 }, { unique: true });

db.images.createIndex({ package: 1, status: 1, queued_at: 1 });  // fila de trabalho e progresso de cada pacote
db.images.createIndex({ package: 1, original_name: 1 });         // import: casar a linha do CSV com a imagem
db.images.createIndex({ equipment: 1, captured_at: -1 });
db.images.createIndex({ camera: 1, captured_at: -1 });
db.images.createIndex({ day_time: 1, captured_at: -1 });
db.images.createIndex({ weather: 1, captured_at: -1 });

db.image_history.createIndex({ image_id: 1, changed_at: -1 });

// Caixas (bounding boxes) do CVAT, uma entrada por tarefa e imagem:
// { task, image: sha1, package, width, height, boxes: [...] }.
// O backend também cria estes índices ao subir (para bancos criados antes dele).
db.createCollection("image_boxes");
db.image_boxes.createIndex({ task: 1, image: 1 }, { unique: true });
db.image_boxes.createIndex({ image: 1 });
db.image_boxes.createIndex({ package: 1 });

// Tarefas de anotação de caixas (ex.: capacete, placa-letras). As caixas de
// image_boxes são por (task, image). O backend também cria este índice ao subir.
db.createCollection("annotation_tasks");
db.annotation_tasks.createIndex({ name: 1 }, { unique: true });

// Perfis de export (os padrões, rf-detr e yolo, ficam no código).
// O backend também cria este índice ao subir.
db.createCollection("export_profiles");
db.export_profiles.createIndex({ name: 1 }, { unique: true });
