import json
from web3 import Web3
from web3 import HTTPProvider
from subprocess import check_output

"""
https://github.com/foundry-rs/foundry

- Init new Project: forge init
- testing: forge test -vvv
"""

RPC_URL = ""
PRIVKEY = ""
SETUP_CONTRACT_ADDR = ""

def get_abi(filename):
    # get abi "solc <filename> --abi"
    abi_str = check_output(['solc', filename, '--abi']).decode().split("Contract JSON ABI")[-1].strip()
    return json.loads(abi_str)

class Account:
    def __init__(self) -> None:
        self.w3 = Web3(HTTPProvider(RPC_URL))
        self.w3.eth.default_account = self.w3.eth.account.from_key(PRIVKEY).address
        self.account_address = self.w3.eth.default_account

    def get_balance(s, addr):
        print("balance:",s.w3.eth.get_balance(addr))


class BaseContract(Account):
    def __init__(self, addr, file, abi=None) -> None:
        super().__init__()
        self.file = file
        self.address = addr
        if abi:
            self.contract = self.w3.eth.contract(addr, abi=abi)
        else:
            self.contract = self.w3.eth.contract(addr, abi=self.get_abi())

    def get_abi(self):
        return get_abi(self.file)


class SetupContract(BaseContract):
    def __init__(self) -> None:
        super().__init__(
            addr=SETUP_CONTRACT_ADDR,
            file="Setup.sol",
        )
    def is_solved(s):
        result = s.contract.functions.isSolved().call()
        print("is solved:", result)

if __name__ == "__main__":
    setup = SetupContract()
    challenge_addr = setup.contract.functions.TARGET().call()
    challenge = ChallengeContract(challenge_addr)
