const fetchData = require('./actual.js');

(async () => {
  try {
    const params = JSON.parse(process.argv[2]);
    console.error(params)
    const data = await fetchData(params.serverURL, params.serverPassword, params.budgetSyncId, params.budgetEncryptionPassword, params.groupName, params.included);

    process.stdout.write(String(JSON.stringify(data) + '\n'))
  } catch (err) {
    console.error('Error: ', err.message || err)
    process.exit(1);
  }
})();
