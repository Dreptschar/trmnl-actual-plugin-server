let api = require('@actual-app/api');
const fs = require('fs');
const path = require("path");

const fetchData = async (serverurl, serverpassword, budgetSyncId, budgetEncPw, groupName, included) => {
    console.error('FETCHING DATA')
    const cacheRootPath = path.resolve('./tmp/cache');
    let folderPath = '';
    let apiInitialised = false;
    try {
        if (typeof budgetSyncId !== 'string' || budgetSyncId.trim() === '') {
            throw new Error('budgetSyncId must be a non-empty string');
        }
        if (budgetEncPw != null && typeof budgetEncPw !== 'string') {
            throw new Error('budgetEncPw must be a string when provided');
        }
        if (typeof groupName !== 'string' || groupName.trim() === '') {
            throw new Error('groupName must be a non-empty string');
        }
        await fs.promises.mkdir(cacheRootPath, { recursive: true });
        folderPath = await fs.promises.mkdtemp(path.join(cacheRootPath, 'run-'));

        await api.init({
            // Budget data will be cached locally here, in subdirectories for each file.
            // This is the URL of your running server
            dataDir: folderPath,
            serverURL: serverurl,
            // This is the password you use to log into the server
            password: serverpassword,
            verbose: true
        });
        apiInitialised = true;
        let mappedCategories = {}
        await api.downloadBudget(
            budgetSyncId,
            budgetEncPw ? { password: budgetEncPw } : undefined
        );
        await api.sync();
        const now = new Date();
        const year = now.getFullYear();
        const month = String(now.getMonth() + 1).padStart(2, '0'); // getMonth() is 0-based
        const formatted = `${year}-${month}`;
        let budget = await api.getBudgetMonth(formatted);
        let categories = budget.categoryGroups.filter(g => g.name === groupName)[0].categories
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
        return { data: mappedCategories };
    } catch (error) {
        console.error("WE CATCHED IT")
        console.error(error)
        return { error: "There was a exception with loading the budget: " + JSON.stringify(serializeErr(error)) }
    } finally {
        try {
            if (apiInitialised) {
                console.error('shuting down')
                await api.shutdown();
            }
        } catch (error) {
            console.error(error)
        }
        try {
            console.error('clearing cache')
            await removeDir(folderPath)
        } catch (error) {
            console.error(error)
        }
    }
}

async function removeDir(dirPath) {
    if (!dirPath) return;
    console.error("Removing cache dir")
    await fs.promises.rm(dirPath, { recursive: true, force: true })
}

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

module.exports = fetchData 
