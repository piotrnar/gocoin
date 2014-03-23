Downloader is a tool designed to quickly download a blockchina from bitcoin network.

You need to start it with the IP of a seed node - some fast hub close to you in the
netowrk is advised. To specify the IP of the seed node use the switch "-s <ip".

The downloader will use the seed node to download all the block headres and then
connect to other peers in order to download the content of the acctual blocks.
As the blocks are being downloaded and parsed the UTXO database is being updated.