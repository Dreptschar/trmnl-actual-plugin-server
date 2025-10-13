const express = require('express');
const fetchData = require('./actual');
const app = express();
const PORT = process.env.PORT || 3000;
app.use(express.json());
app.use(express.urlencoded({ extended: true }));
let locked = false;
// Keep the process alive on late rejections/exceptions
process.on('unhandledRejection', (reason) => {
    console.error('[unhandledRejection]', reason);
});
process.on('uncaughtException', (err) => {
    console.error('[uncaughtException]', err);
    // don't throw here; you want the server to keep running
});
async function withLock(fn) {
    while (locked) await new Promise(r => setTimeout(r, 10));
    locked = true;
    try { return await fn(); }
    finally { locked = false; }
}
app.post('/api/markup', async (req, res, next) => {
    try {
        const body = req.body;
        const data = await withLock(() => fetchData(body.serverURL, body.serverPassword, body.budgetSyncId, body.budgetEncryptionPassword, body.groupName, body.included));
        res.status(200).json(data);
    } catch (err) {
        // Even if something blew up outside fetchData, still 200
        res.status(200).json({ error: JSON.stringify(serializeErr(err)) });
    }
});

function serializeErr(e) {
    if (!e) return null;
    if (e instanceof Error) {
        return {
            name: e.name,
            message: e.message,
            code: e.code,
            stack: e.stack,
        };
    }
    // Non-Error rejections
    return { message: String(e) };
}

app.listen(PORT, () => {
    console.log(`Server listening on port ${PORT}`);
});
