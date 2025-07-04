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

async function fetchData(serverurl, serverpassword, budgetSyncId, budgetEncPw) {
  let api = await require('@actual-app/api');
  const fs = await require('fs');
  const folderPath = '/tmp/cache';
  if (!fs.existsSync(folderPath)) {
    fs.mkdirSync(folderPath, { recursive: true });
    console.log('Folder created:', folderPath);
  }
  await api.init({
    // Budget data will be cached locally here, in subdirectories for each file.
    dataDir: folderPath,
    // This is the URL of your running server
    serverURL: serverurl,
    // This is the password you use to log into the server
    password: serverpassword,
  });
  if(budgetEncPw == null){
  await api.downloadBudget(budgetSyncId);
  }else{
  await api.downloadBudget(budgetSyncId, {
    password: budgetEncPw,
  });
  }
const now = new Date();
const year = now.getFullYear();
const month = String(now.getMonth() + 1).padStart(2, '0'); // getMonth() is 0-based

const formatted = `${year}-${month}`;

  let budget = await api.getBudgetMonth(formatted);
  let categories = budget.categoryGroups[1].categories
  await api.shutdown();

  console.log(budget);
  console.log(categories)

  return categories;
}
