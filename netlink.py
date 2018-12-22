import pprint
import signal
import sys
from enum import Enum

from pyroute2 import IPDB

# http://fos.tech/posts/pyroute2-linux-networking-made-easy/


class NetlinkEvents(Enum):
    # A new neighbor has appeared
    RTM_NEWNEIGH = 'RTM_NEWNEIGH'
    # We're no longer watching a certain neighbor
    RTM_DELNEIGH = 'RTM_DELNEIGH'
    # A new network interface has been created
    RTM_NEWLINK = 'RTM_NEWLINK'
    # A network interface has been deleted
    RTM_DELLINK = 'RTM_DELLINK'
    # An IP address has been added to a network interface
    RTM_NEWADDR = 'RTM_NEWADDR'
    # An IP address has been deleted off of a network interface
    RTM_DELADDR = 'RTM_DELADDR'
    # A route has been added to the routing table
    RTM_NEWROUTE = 'RTM_NEWROUTE'
    # A route has been removed from the routing table
    RTM_DELROUTE = 'RTM_DELROUTE'


ipdb = IPDB()
pp = pprint.PrettyPrinter(indent=3)


def new_event_callback(ipdb, netlink_message, action):
    if True:  # action == NetlinkEvents.RTM_NEWADDR.name:
        print action
        pp.pprint(netlink_message)
        print 100*'-'


if __name__ == "__main__":

    event_callback = ipdb.register_callback(new_event_callback)

    def cleanup(*args):
        ipdb.unregister_callback(event_callback)
        sys.exit()

    signal.signal(signal.SIGINT, cleanup)
    signal.signal(signal.SIGTERM, cleanup)
    signal.pause()
