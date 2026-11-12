// Cria o usuário da aplicação com acesso apenas ao banco sidm.
// Roda só na PRIMEIRA criação do volume, autenticado como root.
db.getSiblingDB("sidm").createUser({
  user: process.env.MONGO_APP_USER,
  pwd: process.env.MONGO_APP_PASSWORD,
  roles: [{ role: "readWrite", db: "sidm" }],
});
