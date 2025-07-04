let api = require('@actual-app/api');

export async function fetchData(serverurl, serverpassword, budgetSyncId, budgetEncPw) {

  await api.init({
    // Budget data will be cached locally here, in subdirectories for each file.
    dataDir: './data/cache',
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
