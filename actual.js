  let api = require('@actual-app/api');
  const fs = require('fs');

const fetchData = async (serverurl, serverpassword, budgetSyncId, budgetEncPw,groupName,excluded) =>{
  const folderPath = '/tmp/cache';
  if (!fs.existsSync(folderPath)) {
    fs.mkdirSync(folderPath, { recursive: true });
  }
  await api.init({
    // Budget data will be cached locally here, in subdirectories for each file.
    // This is the URL of your running server
    dataDir: folderPath,
    serverURL: serverurl,
    // This is the password you use to log into the server
    password: serverpassword,
  });
  let mappedCategories = {}
  try{
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
  let categories = budget.categoryGroups.filter(g => g.name === groupName)[0].categories
  await api.shutdown();
  mappedCategories = categories.filter((x) => !x.hidden).map((c)=> ({ 
    name: c.name,
    budgeted: api.utils.integerToAmount(c.budgeted),
    spent: api.utils.integerToAmount(c.spent),
    balance: api.utils.integerToAmount(c.balance)

  }))
  if(excluded != null){
    const input = excluded;
    const excludedCategories = input.split(",").map(i => i.trim());
    mappedCategories = mappedCategories.filter(c => !excludedCategories.includes(c.name.trim()))
  }
  }catch {
    api.shutdown();
  }

  return mappedCategories;
}

module.exports = fetchData 