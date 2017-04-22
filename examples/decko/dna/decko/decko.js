// Get list cards with chains
expose("listCards", HC.JSON);
function listChains() {return getlink(App.DNA.Hash, "posts");}

// Authorize a new agent_id to participate in this holochain
// agent_id must match the string they use to "hc init" their holochain, and is currently their email by convention
expose("addCard", HC.STRING);
function addMember(x) {
  putmeta(App.DNA.Hash, x, "post")
}

// Initialize by adding agent to holochain
function genesis() {
  //putmeta(App.DNA.Hash, App.Agent.Hash, "post");
  //putmeta(App.DNA.Hash, App.Agent.Hash, "post");
  return true;
}

function validatePut(entry_type,entry,header,sources) {
    return validate(entry_type,entry,header,sources);
}
function validateCommit(entry_type,entry,header,sources) {
    return validate(entry_type,entry,header,sources);
}
// Local validate an entry before committing ???
function validate(entry_type,entry,header,sources) {
    return false;
}

function validateLink(linkingEntryType,baseHash,linkHash,tag,sources){return true}
