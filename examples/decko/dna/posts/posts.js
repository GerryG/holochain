// Get list of posts in a Space
expose("listPosts", HC.JSON);
function listPosts(name) {
  var posts = getlink(name, "posts");
  if( posts instanceof Error ) {
    return []
  } else {
    posts = posts.Entries
    var return_posts = new Array(posts.length);
    for( i=0; i<posts.length; i++) {
      return_posts[i] = JSON.parse(posts[i]["E"]["C"])
      return_posts[i].id = posts[i]["H"]
    }
    return return_posts
  }
}

// Post an event to a card (one that has been newCard(c)'ed already)
expose("postToCard", HC.JSON);
function postToCard(card) {
  card.updateTimestamp = new Date();
  var key = commit("post", card);
  put(key)
  putmeta(card.post, key, "post")
  return key
}

// Create a new card/holochain interface
expose("newCard", HC.JSON);
function newCard(card) {
  card.updateTimestamp = new Date();
  var key = commit("post", card);
  put(key)
  putmeta(card.post, key, "post")
  return key
}

function genesis() {
  return true;
}

function validatePut(entry_type,entry,header,sources) {
    return validate(entry_type,entry,header,sources);
}
function validateCommit(entry_type,entry,header,sources) {
    return validate(entry_type,entry,header,sources);
}

function isValidPost(post) {
    //if !exists(post.name) {
    //	newCard(post)
    //}
    postToCard(post)
    return true
}
// Local validate an entry before committing
function validate(entry_type,entry,header,sources) {
    if( !isValidPost(entry.post) ) {
        debug("post not valid because post "+entry.post.name+" could not be added");
        return false;
    }
    postToCard(post)
}
function validateLink(linkingEntryType,baseHash,linkHash,tag,sources){return true}
