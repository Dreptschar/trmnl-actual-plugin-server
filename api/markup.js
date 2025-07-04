export default function handler(req, res) {
  if (req.method === 'POST') {
    const data = {"text":"Random text 85 from web server","author":"TRMNL","collection":[{"title":"Book title","description":"Some description"},{"title":"Another book","description":"Another description"}]}
    res.status(200).json(JSON.stringify(data));
  } else {
    res.status(405).json({ message: 'Method Not Allowed' });
  }
}