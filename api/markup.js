import { fetchData } from "../actual/actual-service";

export default async function handler(req, res) {
  if (req.method === 'POST') {
    const body = req.body;
    console.log(body)
    const data = await fetchData(body.serverURL, body.serverPassword, body.budgetSyncId, body.budgetEncryptionPassword);
    res.status(200).json(JSON.stringify(data));
  } else {
    res.status(405).json({ message: 'Method Not Allowed' });
  }
}