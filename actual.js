let api = require('@actual-app/api');
const fs = require('fs');
const path = require("path");

const fetchData = async (serverurl, serverpassword, budgetSyncId, budgetEncPw, groupName, included) => {
    const folderPath = './tmp/cache';
    if (!fs.existsSync(folderPath)) {
        fs.mkdirSync(folderPath, {recursive: true});
    }
    await api.init({
        // Budget data will be cached locally here, in subdirectories for each file.
        // This is the URL of your running server
        dataDir: folderPath,
        serverURL: serverurl,
        // This is the password you use to log into the server
        password: serverpassword,
        verbose: true
    });


    let mappedCategories = {}
    try {
        if (budgetEncPw == null) {
            await api.downloadBudget(budgetSyncId);
        } else {
            await api.downloadBudget(budgetSyncId, {
                password: budgetEncPw,
            });
        }
        await api.sync();
        const now = new Date();
        const year = now.getFullYear();
        const month = String(now.getMonth() + 1).padStart(2, '0'); // getMonth() is 0-based

        const formatted = `${year}-${month}`;
        let budget = await api.getBudgetMonth(formatted);
        let categories = budget.categoryGroups.filter(g => g.name === groupName)[0].categories
        await api.shutdown();
        await cleanCache(folderPath)
        mappedCategories = categories.filter((x) => !x.hidden).map((c) => ({
            name: c.name,
            budgeted: api.utils.integerToAmount(c.budgeted),
            spent: api.utils.integerToAmount(c.spent),
            balance: api.utils.integerToAmount(c.balance)

        }))
        if (included != null) {
            const input = included;
            const includedCategories = input.split(",").map(i => i.trim());
            mappedCategories = mappedCategories.filter(c => includedCategories.includes(c.name.trim()))
        }
    } catch(error) {
        await api.shutdown();
        await cleanCache(folderPath)
        return {error: "There was a exception with loading the budget: " + JSON.stringify(error)}
    }

    return mappedCategories;
}

async function cleanCache(dirPath) {
    const entries = await fs.promises.readdir(dirPath)
    console.log(entries)
    for (const entry of entries){
        const fullPath = path.join(dirPath,entry);
        await fs.promises.rm(fullPath, {recursive: true, force: true})
    }
}

module.exports = fetchData 