  let api = require('@actual-app/api');
  const fs = require('fs');

const fetchData = async (serverurl, serverpassword, budgetSyncId, budgetEncPw) =>{
  const folderPath = '/tmp/cache';
  if (!fs.existsSync(folderPath)) {
    fs.mkdirSync(folderPath, { recursive: true });
    console.log('Folder created:', folderPath);
  }
  await api.init({
    // Budget data will be cached locally here, in subdirectories for each file.
    // This is the URL of your running server
    dataDir: folderPath,
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

module.exports = fetchData
