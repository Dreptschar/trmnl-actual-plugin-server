const express = require('express');
const fetchData = require('./actual');
const app = express();
const PORT = process.env.PORT || 3000;

app.use(express.json());

app.post('/api/markup',async (req, res)  => {
    const body = req.body;
    console.log(body)
    const data = await fetchData(body.serverURL, body.serverPassword, body.budgetSyncId, body.budgetEncryptionPassword);
    res.status(200).json(data);
});

app.listen(PORT, () => {
  console.log(`Server listening on port ${PORT}`);
});