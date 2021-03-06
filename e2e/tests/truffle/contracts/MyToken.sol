pragma solidity ^0.4.24;

import "openzeppelin-solidity/contracts/token/ERC721/ERC721Token.sol";

contract MyToken is ERC721Token {
    constructor() ERC721Token("MyToken", "MTC") public {
    }

    function mintToken(uint256 _uid) public {
        _mint(msg.sender, _uid);
    }

    // Workaround for Truffle v4 not handling safeTransferFrom overloads correctly
    function transferToken(address _receiver, uint256 _uid) public {
        safeTransferFrom(msg.sender, _receiver, _uid);
    }
}
